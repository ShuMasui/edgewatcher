package com.edgewatcher.di;

import android.content.Context;
import com.edgewatcher.infrastructure.db.ObservationDatabase;
import dagger.internal.DaggerGenerated;
import dagger.internal.Factory;
import dagger.internal.Preconditions;
import dagger.internal.Provider;
import dagger.internal.QualifierMetadata;
import dagger.internal.ScopeMetadata;
import javax.annotation.processing.Generated;

@ScopeMetadata("javax.inject.Singleton")
@QualifierMetadata("dagger.hilt.android.qualifiers.ApplicationContext")
@DaggerGenerated
@Generated(
    value = "dagger.internal.codegen.ComponentProcessor",
    comments = "https://dagger.dev"
)
@SuppressWarnings({
    "unchecked",
    "rawtypes",
    "KotlinInternal",
    "KotlinInternalInJava",
    "cast",
    "deprecation",
    "nullness:initialization.field.uninitialized"
})
public final class AppModule_DatabaseFactory implements Factory<ObservationDatabase> {
  private final Provider<Context> contextProvider;

  private AppModule_DatabaseFactory(Provider<Context> contextProvider) {
    this.contextProvider = contextProvider;
  }

  @Override
  public ObservationDatabase get() {
    return database(contextProvider.get());
  }

  public static AppModule_DatabaseFactory create(Provider<Context> contextProvider) {
    return new AppModule_DatabaseFactory(contextProvider);
  }

  public static ObservationDatabase database(Context context) {
    return Preconditions.checkNotNullFromProvides(AppModule.INSTANCE.database(context));
  }
}
